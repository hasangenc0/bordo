# ADR-0002: Monorepo + Go Workspace

**Status:** Accepted  
**Date:** 2026-09-26  
**Deciders:** Project founders

## Context

Bordo spans multiple components: control plane, CLI, fleet agent, build layer,
release layer, observe layer, AI agent, web console, Java app frameworks, TS frameworks,
and golden-path templates. We need to decide how to organize the source code.

## Decision

Single git repository (monorepo) with one top-level `go.work` workspace covering all
Go modules. Each major component is a separate Go module in its own directory.
Non-Go components (console, Java SDK, Node SDK) live in their own directories within
the same repo with their own package manager files (`package.json`, `pom.xml`).

```
bordo/
  go.work                  ← covers control-plane, cli, fleet, build, release, observe, agent
  control-plane/go.mod
  cli/go.mod
  fleet/go.mod
  ...
  console/package.json     ← separate; not in go.work
  sdk/java/pom.xml         ← separate
  sdk/node/package.json    ← separate
```

## Alternatives considered

- **Separate repos per component** — better isolation, but makes atomic cross-component
  changes harder, complicates issue tracking, and adds overhead for a small team.
- **Single Go module at root** — simpler initially, but leads to a giant import graph
  with no component boundaries enforced at the module level.
- **Polyrepo with a meta-repo** — too much meta-tooling for the project's current scale.

## Consequences

- Cross-component refactors can be done in a single PR.
- `go work sync` keeps module dependencies consistent.
- GitHub Actions can scope jobs to changed components using path filters.
- Components can still be versioned and released independently by tagging
  `control-plane/v0.1.0` etc. (Go modules support this).
- The repo will grow large; we accept that and manage it with good `.gitignore` and
  sparse-checkout guidance for contributors who only work on one component.
