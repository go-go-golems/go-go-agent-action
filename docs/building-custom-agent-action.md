---
Title: Building and Integrating a Custom Go Review Action
Slug: build-custom-agent-action
Short: Step-by-step guide for cloning the Go agent review template, wiring your own reviewer, and integrating it into GitHub workflows.
Topics:
- github-actions
- code-review
- go
IsTemplate: false
IsTopLevel: false
ShowPerDefault: true
SectionType: GeneralTopic
---

# Building and Integrating a Custom Go Review Action

This tutorial walks you through cloning the Go agent action template, replacing the mock reviewer with your own binary or service, and deploying the action in GitHub workflows. Follow the steps to tailor automated pull-request reviews to your organisation.

## Prerequisites

You should be comfortable with Go modules, Docker container actions, and Git/GitHub basics. Install Go 1.22 or newer and Docker Desktop (or an equivalent engine) locally so you can run unit tests and container builds.

## Step 1 – Clone the template action

Start from the published repository and rename the module to match your GitHub namespace.

```bash
# Fork or copy the template
git clone git@github.com:go-go-golems/go-go-agent-action.git my-agent-action
cd my-agent-action

# Update the module path and imports
perl -pi -e 's#github.com/go-go-golems/go-go-agent-action#github.com/your-org/my-agent-action#g' go.mod cmd/agent-action/main.go
```

Run `GOCACHE=$(pwd)/.cache go test ./...` to ensure the renamed module still builds. This catches typos before you add custom code.

## Step 2 – Implement your reviewer

The action expects a `ReviewResult` JSON from either `tool_mode=http` or `tool_mode=cmd`. Choose the mode that best matches your backend.

### Option A: HTTP service

If you already have a service that generates review feedback, expose a POST endpoint that accepts `PRContext` JSON and emits `ReviewResult`. Configure the action with:

```yaml
with:
  tool_mode: http
  tool_url: https://reviewer.internal/api/review
  tool_token: ${{ secrets.REVIEW_TOKEN }}
  include_patch: true
  output_mode: review+summary
```

### Option B: CLI reviewer

To ship a binary inside the action image, add a new package (for example `cmd/custom-reviewer`) that reads stdin and writes `ReviewResult` to stdout. Wire it in `cmd/agent-action/main.go` so `tool_mode: custom` selects your binary:

```go
case "custom":
    return &action.CommandTool{Command: "/usr/local/bin/custom-reviewer"}, nil
```

Update `Dockerfile` to build and copy the binary into the runtime image. Run `act` to confirm the container invokes your reviewer correctly.

## Step 3 – Configure inputs and triggers

Expose any new knobs in `action.yml`. For example, add `reviewer_config_path` if the reviewer needs a YAML file. Update `internal/action/config.go` to parse the input, and load the file before calling the reviewer. Common patterns include:

- Extending `internal/action/context.go` to add extra metadata (e.g. test results).
- Adjusting `internal/action/triggers.go` to support labels, branches, or comment phrases.
- Adding new `output_mode` combinations in `internal/action/publisher.go` if you want to post to external services.

Document the new inputs in the README so downstream consumers know how to configure them.

## Step 4 – Test locally

Use unit tests and `act` to exercise both pull-request and mention triggers.

```bash
GOCACHE=$(pwd)/.cache go test ./...
~/go/bin/act -W examples/review.yml -P ubuntu-latest=ghcr.io/catthehacker/ubuntu:act-latest
```

Populate `examples/review.yml` with the workflow you expect downstream teams to use. Include `github_token: ${{ secrets.GITHUB_TOKEN }}` so the review can post comments during local simulation.

## Step 5 – Publish the action

1. Commit your changes and push to GitHub.
2. Tag a release (`git tag v1.0.0 && git push origin v1.0.0`).
3. Confirm the tag appears under **Releases** and `action.yml` references the local Dockerfile.

Downstream workflows can now reference `uses: your-org/my-agent-action@v1.0.0`.

## Step 6 – Integrate in a repository

Create a workflow similar to the one `go-go-labs` uses after the merger of PR and mention triggers.

```yaml
name: go-agent-review

on:
  pull_request:
    types: [opened, synchronize, reopened]
  issue_comment:
    types: [created]

permissions:
  contents: read
  pull-requests: write

jobs:
  review:
    if: |
      github.event_name == 'pull_request' ||
      (github.event_name == 'issue_comment' && github.event.issue.pull_request && contains(github.event.comment.body, '@agent'))
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          persist-credentials: false

      - name: Build reviewer
        run: |
          mkdir -p .tooling
          go build -o .tooling/custom-reviewer ./cmd/reviewers/custom-reviewer

      - name: Automated review
        uses: your-org/my-agent-action@v1.0.0
        with:
          tool_mode: cmd
          tool_cmd: ./.tooling/custom-reviewer
          include_patch: true
          output_mode: review+summary
          github_token: ${{ secrets.GITHUB_TOKEN }}
```

Developers receive automated reviews every time they update a PR or explicitly call the reviewer with `@agent`. Because the reviewer binary lives in the repository under test, you can evolve scripts and configuration alongside the application code.

## Step 7 – Extend and iterate

As your review needs grow, consider:

- Allowing reviewers to run optional suites by passing structured commands through the action inputs.
- Aggregating results from multiple tools (lint, security scan, QA scripts) before posting a single set of comments.
- Publishing a Checks API summary or custom artifacts from `internal/action/publisher.go` for richer CI dashboards.

Each enhancement should come with updated documentation, examples, and ideally automated tests that you can run with `act` or unit suites.

## Troubleshooting tips

- **401 errors when creating reviews** – Ensure `github_token` (or a PAT) is passed in workflow inputs. The default `mock` mode cannot post without it.
- **422 “diff hunk” errors** – Avoid inline comments when running from `issue_comment` events unless your reviewer knows the diff context. Use issue comments or fall back to the pull-request trigger.
- **Docker build failures** – If BuildKit flags cache mounts, remove the `RUN --mount` directives from the Dockerfile, or enable BuildKit explicitly.

With these steps you can deliver a tailored review experience while reusing the Go agent action’s proven GitHub integration.
