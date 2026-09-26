# Contributing a Golden-Path Template

Templates live in `templates/<name>/`. Follow this guide to add a new one.

## Template directory layout

```
templates/my-template/
  README.md         # required — describes what this template produces
  template.yaml     # required — Bordo template manifest
  Dockerfile        # required for buildable templates
  src/              # source code with {{.Var}} placeholders
  ...
```

## template.yaml

```yaml
# template.yaml
name: my-template
description: One-line description
language: java           # java | typescript | go | other
runtime: web-service     # web-service | worker | kafka | job | frontend | bff | library
variables:
  - name: ProjectName
    prompt: "Project name (kebab-case, e.g. my-service)"
    default: ""
    required: true
  - name: GroupId
    prompt: "Maven group ID (e.g. com.example)"
    default: "com.example"
    required: false
build:
  dockerfile: Dockerfile  # relative to template root
  context: .
health:
  path: /actuator/health  # HTTP path Bordo polls after deploy
  port: 8080
observability:
  metrics: true           # bordo injects OTel sidecar if true
  logs: true
```

## Variable placeholders

Use Go template syntax in any text file:

```
// Package {{.ProjectName}} is the entry point.
package {{.ProjectName | lower | replace "-" "_"}}
```

Available variables:
- `{{.ProjectName}}` — from the `name` variable
- `{{.GroupId}}` — from the `groupId` variable (Java)
- `{{.BordoVersion}}` — the Bordo version that created the project
- `{{.CreatedAt}}` — ISO8601 timestamp

## Testing your template

```bash
# Expand the template locally without registering it
bordo template expand templates/my-template \
  --var ProjectName=test-svc \
  --output /tmp/test-svc

# Build the expanded project
cd /tmp/test-svc && docker build .
```

## PR checklist

- [ ] `template.yaml` passes schema validation (`bordo template validate templates/my-template`)
- [ ] Template expands without errors with test vars
- [ ] Expanded project builds (Dockerfile works)
- [ ] `README.md` describes: what it produces, variables, build output, and any config
- [ ] Health check path is correct and responds 200 after startup
- [ ] Tracker issue `BRD-006` (or a new issue) references this PR
