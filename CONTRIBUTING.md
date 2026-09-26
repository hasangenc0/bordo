# Contributing to Bordo

Thank you for contributing! Bordo is organized as a filesystem kanban board; all work
is tracked in `tracker/`. Read this and `tracker/README.md` before opening a PR.

## Process

1. **Find or create an issue** in `tracker/backlog/` or `tracker/todo/`.
2. **Move it to `in-progress/`** and set `assignee:` to your GitHub handle.
3. **Work on it** in a branch named `brd-<id>-<short-slug>`.
4. **PR targeting `main`** — title: `BRD-<id>: <title>`. CI must pass.
5. **Review**: move issue to `review/`, reference the PR in the notes log.
6. **Merge & done**: move issue to `done/`, set `updated:`.

## Code standards

- **Go**: `gofmt` + `goimports`. Run `make fmt` before committing.
- **Java**: Google Java Format (formatter config in `sdk/java/`).
- **TypeScript**: `prettier` (config at `console/.prettierrc`).
- **Commits**: present tense imperative — `Add project registry`, not `Added`.
- **Secrets**: never commit credentials, tokens, or keys. Use `.env.example` for shapes.

## Adding a golden-path template

Templates live in `templates/<name>/`. Each must include:
- `README.md` — what this template produces, how to use it, config options
- `template.yaml` — Bordo template manifest (schema in `api/openapi/`)
- Source files (with `{{.ProjectName}}` etc. placeholders where appropriate)

See `docs/guides/contributing-a-template.md` for the full contract.

## Running locally

```bash
# Build everything
make build

# Run the control plane (listens :7400 gRPC, :7401 HTTP)
./bin/bordod serve --dev

# Use the CLI against it
./bin/bordo --server localhost:7400 project list
```

## Issue IDs

Do not reuse IDs. Check `tracker/` for the highest existing `BRD-<id>` and increment.
See `tracker/ISSUE_TEMPLATE.md` for the canonical file format.

## License

By contributing you agree your contributions are licensed under Apache 2.0.
