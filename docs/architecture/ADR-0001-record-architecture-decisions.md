# ADR-0001: Record Architecture Decisions

**Status:** Accepted  
**Date:** 2026-09-26  
**Deciders:** Project founders

## Context

As an open-source project with parallel workstreams, we need a lightweight way to
capture significant architectural decisions — including the context and alternatives
considered — so that future contributors can understand why the codebase is shaped
the way it is without having to reconstruct the reasoning.

## Decision

We will use Architecture Decision Records (ADRs), stored as Markdown files in
`docs/architecture/`, named `ADR-NNNN-<short-title>.md`.

Format:
- **Status** — Proposed / Accepted / Deprecated / Superseded-by ADR-XXXX
- **Date**
- **Deciders**
- **Context** — the problem
- **Decision** — what we chose
- **Alternatives considered**
- **Consequences**

ADRs are immutable once accepted. To change a decision, create a new ADR that supersedes it.

## Alternatives considered

- Wiki pages — too easy to silently edit, no history
- GitHub Issues — good for discussion, poor for archival reference
- Inline comments in code — not discoverable enough for cross-cutting decisions

## Consequences

- Contributors have a searchable record of past decisions.
- New ADRs require a short PR — the review process adds intentionality.
- The number grows over time; that is intentional (it's a log, not a clean slate).
